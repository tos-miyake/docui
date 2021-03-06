package panel

import (
	"fmt"
	"os"
	"strings"
	"time"

	docker "github.com/fsouza/go-dockerclient"
	"github.com/jroimartin/gocui"
	"github.com/skanehira/docui/common"
)

type ImageList struct {
	*Gui
	name string
	Position
	Images         []*Image
	Data           map[string]interface{}
	ClosePanelName string
	Items          Items
	selectedImage  *Image
	filter         string
}

type Image struct {
	ID      string `tag:"ID" len:"min:0.1 max:0.2"`
	Repo    string `tag:"REPOSITORY" len:"min:0.1 max:0.3"`
	Tag     string `tag:"TAG" len:"min:0.1 max:0.1"`
	Created string `tag:"CREATED" len:"min:0.1 max:0.2"`
	Size    string `tag:"SIZE" len:"min:0.1 max:0.2"`
}

func NewImageList(gui *Gui, name string, x, y, w, h int) *ImageList {
	i := &ImageList{
		Gui:      gui,
		name:     name,
		Position: Position{x, y, w, h},
		Data:     make(map[string]interface{}),
		Items:    Items{},
	}

	return i
}

func (i *ImageList) Name() string {
	return i.name
}

func (i *ImageList) Edit(v *gocui.View, key gocui.Key, ch rune, mod gocui.Modifier) {
	switch {
	case ch != 0 && mod == 0:
		v.EditWrite(ch)
	case key == gocui.KeySpace:
		v.EditWrite(' ')
	case key == gocui.KeyBackspace || key == gocui.KeyBackspace2:
		v.EditDelete(true)
	case key == gocui.KeyArrowLeft:
		v.MoveCursor(-1, 0, false)
		return
	case key == gocui.KeyArrowRight:
		v.MoveCursor(+1, 0, false)
		return
	}

	i.filter = ReadLine(v, nil)

	if v, err := i.View(i.name); err == nil {
		i.GetImageList(v)
	}
}

func (i *ImageList) SetView(g *gocui.Gui) error {
	// set header panel
	if v, err := g.SetView(ImageListHeaderPanel, i.x, i.y, i.w, i.h); err != nil {
		if err != gocui.ErrUnknownView {
			panic(err)
		}

		v.Wrap = true
		v.Frame = true
		v.Title = v.Name()
		v.FgColor = gocui.AttrBold | gocui.ColorWhite
		common.OutputFormatedHeader(v, &Image{})
	}

	// set scroll panel
	v, err := g.SetView(i.name, i.x, i.y+1, i.w, i.h)
	if err != nil {
		if err != gocui.ErrUnknownView {
			return err
		}
		v.Frame = false
		v.Wrap = true
		v.FgColor = gocui.ColorCyan
		v.SelBgColor = gocui.ColorWhite
		v.SelFgColor = gocui.ColorBlack | gocui.AttrBold
		v.SetOrigin(0, 0)
		v.SetCursor(0, 0)

		i.GetImageList(v)
	}

	i.SetKeyBinding()

	//  monitoring container status interval 5s
	go func() {
		for {
			i.Update(func(g *gocui.Gui) error {
				i.Refresh(g, v)
				return nil
			})
			time.Sleep(5 * time.Second)
		}
	}()

	return nil
}

func (i *ImageList) Refresh(g *gocui.Gui, v *gocui.View) error {
	i.Update(func(g *gocui.Gui) error {
		v, err := i.View(i.name)
		if err != nil {
			panic(err)
		}
		i.GetImageList(v)
		return nil
	})

	return nil
}

func (i *ImageList) SetKeyBinding() {
	i.SetKeyBindingToPanel(i.name)

	if err := i.SetKeybinding(i.name, 'j', gocui.ModNone, CursorDown); err != nil {
		panic(err)
	}
	if err := i.SetKeybinding(i.name, 'k', gocui.ModNone, CursorUp); err != nil {
		panic(err)
	}
	if err := i.SetKeybinding(i.name, gocui.KeyEnter, gocui.ModNone, i.DetailImage); err != nil {
		panic(err)
	}
	if err := i.SetKeybinding(i.name, 'o', gocui.ModNone, i.DetailImage); err != nil {
		panic(err)
	}
	if err := i.SetKeybinding(i.name, 'c', gocui.ModNone, i.CreateContainerPanel); err != nil {
		panic(err)
	}
	if err := i.SetKeybinding(i.name, 'p', gocui.ModNone, i.PullImagePanel); err != nil {
		panic(err)
	}
	if err := i.SetKeybinding(i.name, 'd', gocui.ModNone, i.RemoveImage); err != nil {
		panic(err)
	}
	if err := i.SetKeybinding(i.name, gocui.KeyCtrlD, gocui.ModNone, i.RemoveDanglingImages); err != nil {
		panic(err)
	}
	if err := i.SetKeybinding(i.name, 's', gocui.ModNone, i.SaveImagePanel); err != nil {
		panic(err)
	}
	if err := i.SetKeybinding(i.name, 'i', gocui.ModNone, i.ImportImagePanel); err != nil {
		panic(err)
	}
	if err := i.SetKeybinding(i.name, gocui.KeyCtrlL, gocui.ModNone, i.LoadImagePanel); err != nil {
		panic(err)
	}
	if err := i.SetKeybinding(i.name, gocui.KeyCtrlS, gocui.ModNone, i.SearchImagePanel); err != nil {
		panic(err)
	}
	if err := i.SetKeybinding(i.name, gocui.KeyCtrlR, gocui.ModNone, i.Refresh); err != nil {
		panic(err)
	}
	if err := i.SetKeybinding(i.name, 'f', gocui.ModNone, i.Filter); err != nil {
		panic(err)
	}
}

func (i *ImageList) selected() (*Image, error) {
	v, _ := i.View(i.name)
	_, cy := v.Cursor()
	_, oy := v.Origin()

	index := oy + cy
	length := len(i.Images)

	if index >= length {
		return nil, common.NoImage
	}

	return i.Images[index], nil
}

func (i *ImageList) CreateContainerPanel(g *gocui.Gui, v *gocui.View) error {
	i.NextPanel = i.name

	name, err := i.GetImageName()
	if err != nil {
		i.ErrMessage(err.Error(), i.NextPanel)
		return nil
	}

	i.Data = map[string]interface{}{
		"Image": name,
	}

	maxX, maxY := i.Size()
	x := maxX / 8
	y := maxY / 8
	w := maxX - x
	h := maxY - y

	i.ClosePanelName = CreateContainerPanel
	i.Items = i.NewCreateContainerItems(x, y, w, h)

	handlers := Handlers{
		gocui.KeyEnter: i.CreateContainer,
	}

	NewInput(i.Gui, CreateContainerPanel, x, y, w, h, i.Items, i.Data, handlers)
	return nil
}

func (i *ImageList) CreateContainer(g *gocui.Gui, v *gocui.View) error {
	data, err := i.GetItemsToMap(i.Items)
	if err != nil {
		i.ClosePanel(g, v)
		i.ErrMessage(err.Error(), i.NextPanel)
		return nil
	}

	options, err := i.Docker.NewContainerOptions(data)

	if err != nil {
		i.ClosePanel(g, v)
		i.ErrMessage(err.Error(), i.NextPanel)
		return nil
	}

	g.Update(func(g *gocui.Gui) error {
		i.ClosePanel(g, v)
		i.StateMessage("container creating...")

		g.Update(func(g *gocui.Gui) error {
			defer i.CloseStateMessage()

			if err := i.Docker.CreateContainerWithOptions(options); err != nil {
				i.ErrMessage(err.Error(), i.NextPanel)
				return nil
			}

			i.Panels[ContainerListPanel].Refresh(g, v)
			i.SwitchPanel(i.NextPanel)

			return nil
		})

		return nil
	})

	return nil
}

func (i *ImageList) PullImagePanel(g *gocui.Gui, v *gocui.View) error {
	maxX, maxY := i.Size()
	x := maxX / 8
	y := maxY / 3
	w := maxX - x
	h := y + 4

	i.NextPanel = i.name
	i.ClosePanelName = PullImagePanel
	i.Items = i.NewPullImageItems(x, y, w, h)

	handlers := Handlers{
		gocui.KeyEnter: i.PullImage,
	}

	NewInput(i.Gui, PullImagePanel, x, y, w, h, i.Items, i.Data, handlers)
	return nil
}

func (i *ImageList) PullImage(g *gocui.Gui, v *gocui.View) error {

	item := strings.SplitN(ReadLine(v, nil), ":", 2)

	if len(item) == 0 {
		return nil
	}

	name := item[0]
	var tag string

	if len(item) == 1 {
		tag = "latest"
	} else {
		tag = item[1]
	}

	g.Update(func(g *gocui.Gui) error {
		i.ClosePanel(g, v)
		i.StateMessage("image pulling...")

		g.Update(func(g *gocui.Gui) error {
			defer i.CloseStateMessage()

			options := docker.PullImageOptions{
				Repository: name,
				Tag:        tag,
			}

			if err := i.Docker.PullImageWithOptions(options); err != nil {
				i.ErrMessage(err.Error(), i.NextPanel)
				return nil
			}

			i.Refresh(g, v)
			i.SwitchPanel(i.NextPanel)

			return nil

		})

		return nil
	})

	return nil
}

func (i *ImageList) DetailImage(g *gocui.Gui, v *gocui.View) error {
	i.NextPanel = i.name

	image, err := i.selected()
	if err != nil {
		i.ErrMessage(err.Error(), i.NextPanel)
		return nil
	}

	img, err := i.Docker.InspectImage(image.ID)
	if err != nil {
		i.ErrMessage(err.Error(), i.NextPanel)
		return nil
	}

	i.PopupDetailPanel(g, v)

	v, err = g.View(DetailPanel)
	if err != nil {
		panic(err)
	}

	v.Clear()
	v.SetOrigin(0, 0)
	v.SetCursor(0, 0)
	fmt.Fprint(v, common.StructToJson(img))

	return nil
}

func (i *ImageList) SaveImagePanel(g *gocui.Gui, v *gocui.View) error {
	i.NextPanel = i.name

	name, err := i.GetImageName()
	if err != nil {
		i.ErrMessage(err.Error(), i.NextPanel)
		return nil
	}

	maxX, maxY := i.Size()
	x := maxX / 8
	y := maxY / 3
	w := maxX - x
	h := y + 4

	i.ClosePanelName = SaveImagePanel
	i.Items = i.NewSaveImageItems(x, y, w, h)

	i.Data = map[string]interface{}{
		"ID": name,
	}

	handlers := Handlers{
		gocui.KeyEnter: i.SaveImage,
	}

	NewInput(i.Gui, SaveImagePanel, x, y, w, h, i.Items, i.Data, handlers)
	return nil
}

func (i *ImageList) SaveImage(g *gocui.Gui, v *gocui.View) error {
	path := ReadLine(v, nil)

	if path == "" {
		return nil
	}

	g.Update(func(g *gocui.Gui) error {
		i.ClosePanel(g, v)
		i.StateMessage("image saving....")

		g.Update(func(g *gocui.Gui) error {
			defer i.CloseStateMessage()

			file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0666)
			if err != nil {
				i.ErrMessage(err.Error(), i.NextPanel)
				return nil
			}
			defer file.Close()

			options := docker.ExportImageOptions{
				Name:         i.Data["ID"].(string),
				OutputStream: file,
			}

			if err := i.Docker.SaveImageWithOptions(options); err != nil {
				i.ErrMessage(err.Error(), i.NextPanel)
				return nil
			}

			i.SwitchPanel(i.NextPanel)

			return nil
		})

		return nil
	})

	return nil
}

func (i *ImageList) ImportImagePanel(g *gocui.Gui, v *gocui.View) error {

	maxX, maxY := i.Size()
	x := maxX / 8
	y := maxY / 3
	w := maxX - x
	h := maxY - y

	i.NextPanel = i.name
	i.ClosePanelName = ImportImagePanel
	i.Items = i.NewImportImageItems(x, y, w, h)

	handlers := Handlers{
		gocui.KeyEnter: i.ImportImage,
	}

	NewInput(i.Gui, ImportImagePanel, x, y, w, h, i.Items, i.Data, handlers)
	return nil
}

func (i *ImageList) ImportImage(g *gocui.Gui, v *gocui.View) error {
	data, err := i.GetItemsToMap(i.Items)
	if err != nil {
		i.ClosePanel(g, v)
		i.ErrMessage(err.Error(), i.NextPanel)
		return nil
	}

	options := docker.ImportImageOptions{
		Repository: data["Repository"],
		Source:     data["Path"],
		Tag:        data["Tag"],
	}

	g.Update(func(g *gocui.Gui) error {
		i.ClosePanel(g, v)
		i.StateMessage("image importing....")

		g.Update(func(g *gocui.Gui) error {
			defer i.CloseStateMessage()

			if err := i.Docker.ImportImageWithOptions(options); err != nil {
				i.ErrMessage(err.Error(), i.NextPanel)
				return nil
			}

			i.Refresh(g, v)
			i.SwitchPanel(i.NextPanel)

			return nil
		})

		return nil
	})

	return nil
}

func (i *ImageList) LoadImagePanel(g *gocui.Gui, v *gocui.View) error {
	i.NextPanel = i.name

	maxX, maxY := i.Size()
	x := maxX / 8
	y := maxY / 3
	w := maxX - x
	h := y + 4

	i.NextPanel = i.name
	i.ClosePanelName = LoadImagePanel
	i.Items = i.NewLoadImageItems(x, y, w, h)

	handlers := Handlers{
		gocui.KeyEnter: i.LoadImage,
	}

	NewInput(i.Gui, LoadImagePanel, x, y, w, h, i.Items, i.Data, handlers)
	return nil
}

func (i *ImageList) LoadImage(g *gocui.Gui, v *gocui.View) error {
	path := ReadLine(v, nil)
	if path == "" {
		return nil
	}

	g.Update(func(g *gocui.Gui) error {
		i.ClosePanel(g, v)
		i.StateMessage("image loading....")

		g.Update(func(g *gocui.Gui) error {

			defer i.CloseStateMessage()
			if err := i.Docker.LoadImageWithPath(path); err != nil {
				i.ErrMessage(err.Error(), i.NextPanel)
				return nil
			}

			i.Refresh(g, v)
			i.SwitchPanel(i.NextPanel)

			return nil
		})

		return nil
	})

	return nil
}

func (i *ImageList) SearchImagePanel(g *gocui.Gui, v *gocui.View) error {
	i.NextPanel = g.CurrentView().Name()

	maxX, maxY := g.Size()
	x := maxX / 8
	y := maxY / 4
	w := maxX - x
	h := y + 2

	NewSearchImage(i.Gui, SearchImagePanel, Position{x, y, w, h})
	return nil
}

func (i *ImageList) GetImageList(v *gocui.View) {
	v.Clear()
	i.Images = make([]*Image, 0)

	for _, image := range i.Docker.Images(docker.ListImagesOptions{}) {
		for _, repoTag := range image.RepoTags {
			repo, tag := ParseRepoTag(repoTag)

			if i.filter != "" {
				name := fmt.Sprintf("%s:%s", repo, tag)
				if strings.Index(strings.ToLower(name), strings.ToLower(i.filter)) == -1 {
					continue
				}
			}

			id := image.ID[7:19]
			created := ParseDateToString(image.Created)
			size := ParseSizeToString(image.Size)

			image := &Image{
				ID:      id,
				Repo:    repo,
				Tag:     tag,
				Created: created,
				Size:    size,
			}

			i.Images = append(i.Images, image)

			common.OutputFormatedLine(v, image)
		}
	}
}

func (i *ImageList) GetImageName() (string, error) {
	image, err := i.selected()
	if err != nil {
		return "", err
	}

	var name string
	if image.Repo == "<none>" || image.Tag == "<none>" {
		name = image.ID
	} else {
		name = fmt.Sprintf("%s:%s", image.Repo, image.Tag)
	}

	return name, nil
}

func (i *ImageList) RemoveImage(g *gocui.Gui, v *gocui.View) error {
	i.NextPanel = i.name

	name, err := i.GetImageName()
	if err != nil {
		i.ErrMessage(err.Error(), i.NextPanel)
		return nil
	}

	i.ConfirmMessage("Are you sure you want to remove this image? (y/n)", func(g *gocui.Gui, v *gocui.View) error {
		defer i.Refresh(g, v)
		defer i.CloseConfirmMessage(g, v)

		if err := i.Docker.RemoveImageWithName(name); err != nil {
			i.ErrMessage(err.Error(), i.NextPanel)
			return nil
		}

		return nil
	})

	return nil
}

func (i *ImageList) RemoveDanglingImages(g *gocui.Gui, v *gocui.View) error {
	i.NextPanel = i.name

	if len(i.Images) == 0 {
		i.ErrMessage(common.NoImage.Error(), i.NextPanel)
		return nil
	}

	i.ConfirmMessage("Are you sure you want to remove dangling images? (y/n)", func(g *gocui.Gui, v *gocui.View) error {
		defer i.Refresh(g, v)
		defer i.CloseConfirmMessage(g, v)

		if err := i.Docker.RemoveDanglingImages(); err != nil {
			i.ErrMessage(err.Error(), i.NextPanel)
			return nil
		}

		return nil
	})
	return nil
}

func (i *ImageList) Filter(g *gocui.Gui, lv *gocui.View) error {
	i.NextPanel = i.name

	isReset := false
	closePanel := func(g *gocui.Gui, v *gocui.View) error {
		if isReset {
			i.filter = ""
		} else {
			lv.SetCursor(0, 0)
			i.filter = ReadLine(v, nil)
		}
		if v, err := i.View(i.name); err == nil {
			i.GetImageList(v)
		}

		if err := g.DeleteView(v.Name()); err != nil {
			panic(err)
		}

		g.DeleteKeybindings(v.Name())
		i.SwitchPanel(i.name)
		return nil
	}

	reset := func(g *gocui.Gui, v *gocui.View) error {
		isReset = true
		return closePanel(g, v)
	}

	if err := i.NewFilterPanel(i, reset, closePanel); err != nil {
		panic(err)
	}

	return nil
}

func (i *ImageList) ClosePanel(g *gocui.Gui, v *gocui.View) error {
	return i.Panels[i.ClosePanelName].(*Input).ClosePanel(g, v)
}

func (i *ImageList) NewSaveImageItems(ix, iy, iw, ih int) Items {
	names := []string{
		"Path",
	}

	return NewItems(names, ix, iy, iw, ih, 6)
}

func (i *ImageList) NewImportImageItems(ix, iy, iw, ih int) Items {
	names := []string{
		"Repository",
		"Path",
		"Tag",
	}

	return NewItems(names, ix, iy, iw, ih, 12)
}

func (i *ImageList) NewLoadImageItems(ix, iy, iw, ih int) Items {
	names := []string{
		"Path",
	}

	return NewItems(names, ix, iy, iw, ih, 6)
}

func (i *ImageList) NewPullImageItems(ix, iy, iw, ih int) Items {
	names := []string{
		"Name",
	}

	return NewItems(names, ix, iy, iw, ih, 6)
}

func (i *ImageList) NewCreateContainerItems(ix, iy, iw, ih int) Items {
	names := []string{
		"Name",
		"HostPort",
		"Port",
		"HostVolume",
		"Volume",
		"Image",
		"Attach",
		"Env",
		"Cmd",
	}

	return NewItems(names, ix, iy, iw, ih, 12)
}
